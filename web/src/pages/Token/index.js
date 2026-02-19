import React from 'react';
import { Container } from 'semantic-ui-react';
import AggTokensTable from '../../components/AggTokensTable';
import Header from '../../components/Header';
import Footer from '../../components/Footer';

const Token = () => (
    <>
        <Header />
        <Container>
            <AggTokensTable />
        </Container>
        <Footer />
    </>
);

export default Token;
