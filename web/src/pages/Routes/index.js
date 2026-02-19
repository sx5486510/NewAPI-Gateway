import React from 'react';
import { Container } from 'semantic-ui-react';
import ModelRoutesTable from '../../components/ModelRoutesTable';
import Header from '../../components/Header';
import Footer from '../../components/Footer';

const Routes = () => (
    <>
        <Header />
        <Container>
            <ModelRoutesTable />
        </Container>
        <Footer />
    </>
);

export default Routes;
